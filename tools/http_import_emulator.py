#! /usr/bin/env python3

# Simulate HTTP import endpoint: accept and display requests.

import argparse
import gzip
import http.client
import http.server
import re
import socketserver
import zlib
from http import HTTPStatus
from typing import BinaryIO, Callable, Optional

DEFAULT_PORT = 8080


class Decoder:
    pass


class DecodeGzip(Decoder):
    def __init__(self, fh: BinaryIO):
        self._fh = gzip.open(fh, "rb")

    def read(self, n: Optional[int] = None) -> bytes:
        return self._fh.read(n)


class DecodeDeflate(Decoder):
    def __init__(self, fh: BinaryIO):
        self._fh = fh
        self._b = None

    def read(self, n: Optional[int] = None) -> bytes:
        if self._b is None:
            self._b = zlib.decompress(self._fh.read())
            self._n_read = 0
            self._content_length = len(self._b)
        if n is None:
            to_read = self._content_length - self._n_read
        else:
            to_read = min(n, self._content_length - self._n_read)
        b = b""
        if to_read > 0:
            if self._n_read == 0 and to_read == self._content_length:
                b = self._b
            else:
                b = self._b[self._n_read : self._n_read + to_read + 1]
            self._n_read += to_read
        return b


SUPPORTED_ENCODING = {
    "gzip": DecodeGzip,
    "x-gzip": DecodeGzip,
    "deflate": DecodeDeflate,
}


class BodyError(Exception):
    pass


class BodyReader:
    def __init__(self, fh: BinaryIO, content_length: int = 0):
        if content_length <= 0:
            raise BodyError(f"{content_length}: Invalid Content-Length")
        self._fh = fh
        self._content_length = content_length
        self._n_read = 0

    def read(self, n: Optional[int] = None) -> bytes:
        if n is None:
            to_read = self._content_length - self._n_read
        else:
            to_read = min(n, self._content_length - self._n_read)
        if to_read > 0:
            b = self._fh.read(to_read)
            self._n_read += len(b)
        else:
            b = b""
        return b


class ChunkReader:
    def __init__(self, fh: BinaryIO, fmt_logger: Optional[Callable] = None):
        self._fh = fh
        self._fmt_logger = fmt_logger
        self._br = None
        self._at_eof = False
        self._locate_next_chunk()

    def _locate_next_chunk(self):
        chunk_sz_ext = b""
        # Read until '\r':
        while True:
            b = self._fh.read(1)
            if len(b) == 0:
                return
            if b == b"\r":
                break
            chunk_sz_ext += b
        # Expect '\n':
        b = self._fh.read(1)
        if b != b"\n":
            return
        # Parse chunk_sz_ext:
        try:
            sz_ext = re.split(r"\s*;\s*", str(chunk_sz_ext, "utf-8"))
            chunk_sz = int(sz_ext[0], base=16)
            chunk_ext = [re.split(r"\s*=\s*", e.strip()) for e in sz_ext[1:]]
        except (ValueError, TypeError, IndexError, UnicodeDecodeError):
            raise BodyError("Cannot process chunk header")
        if self._fmt_logger is not None:
            self._fmt_logger(
                "Chunk size: %d, extensions: %s",
                chunk_sz,
                repr(chunk_ext),
            )
        if chunk_sz > 0:
            self._br = BodyReader(self._fh, chunk_sz)
        else:
            self._verify_end_of_chunk()

    def _verify_end_of_chunk(self):
        if not self._at_eof:
            if self._fh.read(2) != b"\r\n":
                raise BodyError("Cannot locate chunk end")
            self._at_eof = True

    def read(self, n: Optional[int] = None) -> bytes:
        if self._at_eof:
            return b""
        b = self._br.read(n)
        if len(b) == 0:
            self._verify_end_of_chunk()
        return b

    @property
    def at_eof(self):
        return self._at_eof


class ChunkedBodyReader:
    def __init__(
        self,
        fh: BinaryIO,
        decoder_class: Optional[Decoder] = None,
        fmt_logger: Optional[Callable] = None,
    ):
        self._fh = fh
        self._decoder_class = decoder_class
        self._fmt_logger = fmt_logger
        self._cr = None
        self._last_chunk = False

    def read(self, n: Optional[int] = None) -> bytes:
        b = b""
        to_read = n
        while (to_read is None or to_read > 0) and not self._last_chunk:
            if self._cr is None:
                cr = ChunkReader(self._fh, fmt_logger=self._fmt_logger)
                if cr.at_eof:
                    cr = None
                    self._last_chunk = True
                    break
                elif self._decoder_class is not None:
                    cr = self._decoder_class(cr)
                self._cr = cr
            chunk_b = self._cr.read(to_read)
            chunk_n = len(chunk_b)
            if chunk_n > 0:
                b += chunk_b
                if to_read is not None:
                    to_read -= chunk_n
            else:
                self._cr = None
        return b


class HTTPRequestHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.log_message("%s\n%s", self.requestline, self.headers)

        transfer_encodings = set(
            re.split(r"\s*,\s*", self.headers.get("Transfer-Encoding", "").strip())
        )
        transfer_decoder_class = None
        is_chunked = False
        for encoding in transfer_encodings:
            if encoding == "":
                continue
            if encoding == "chunked":
                is_chunked = True
                continue
            transfer_decoder_class = SUPPORTED_ENCODING.get(encoding)
            if transfer_decoder_class is None:
                self.send_error(
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE,
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE.description,
                    explain=f"{encoding}: unsupported Transfer-Encoding",
                )
                return
        if is_chunked:
            body_reader = ChunkedBodyReader(
                self.rfile, transfer_decoder_class, self.log_message
            )
        else:
            try:
                content_length = int(self.headers["Content-Length"])
                body_reader = BodyReader(self.rfile, content_length)
            except (ValueError, TypeError, KeyError, BodyError):
                self.send_error(
                    HTTPStatus.LENGTH_REQUIRED,
                    HTTPStatus.LENGTH_REQUIRED.description,
                    explain="Missing/invalid Content-Length",
                )
                return
            if transfer_decoder_class is not None:
                body_reader = transfer_decoder_class(body_reader)

        content_encodings = re.split(
            r"\s*,\s*", self.headers.get("Content-Encoding", "").strip()
        )
        content_decoder_class = None
        for encoding in content_encodings:
            if encoding == "":
                pass
            content_decoder_class = SUPPORTED_ENCODING.get(encoding)
            if content_decoder_class is None:
                self.send_error(
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE,
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE.description,
                    explain=f"{encoding}: unsupported Content-Encoding",
                )
                return
        if content_decoder_class is not None:
            body_reader = content_decoder_class(body_reader)

        expect_match = re.match("\s*(\d+)", self.headers.get("Expect", ""))
        if expect_match is not None:
            self.send_response(HTTPStatus(int(expect_match.groups()[0])))
            self.end_headers()

        try:
            body = body_reader.read()
        except (BodyError):
            self.send_error(
                HTTPStatus.UNPROCESSABLE_ENTITY,
                HTTPStatus.UNPROCESSABLE_ENTITY.description,
            )
            return
        self.log_message("Body: %d bytes after decoding", len(body))

        if self.server._context.get("show_body"):
            content_type = (
                self.headers.get_content_type() or self.headers.get_default_type()
            )
            content_main_type, content_sub_type = content_type.split("/")
            if content_main_type == "text":
                body = str(body, "utf-8")
            self.log_message("\n%s\n", body)

        self.send_response(HTTPStatus.OK)
        self.end_headers()

    def do_GET(self):
        self.log_message(f""""{self.requestline}"\n{self.headers}""")
        self.send_response(HTTPStatus.OK)
        self.end_headers()

    do_HEAD = do_GET
    do_PUT = do_POST


class HttpServerWithContext(socketserver.ThreadingTCPServer):
    def __init__(self, address_port, handler, **context):
        super().__init__(address_port, handler)
        self._context = context


def run(
    address="",
    port=DEFAULT_PORT,
    **context,
):
    with HttpServerWithContext((address, port), HTTPRequestHandler, **context) as httpd:
        print(f"Accepting connections to {'*' if not address else address}:{port}")
        httpd.serve_forever()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-p",
        "--port",
        type=int,
        default=DEFAULT_PORT,
        help="""
            Port to listen to for connections, default: %(default)d
        """,
    )
    parser.add_argument(
        "-b",
        "--bind-address",
        default="",
        help="""
            Bind to a specific address, default: %(default)s
        """,
    )
    parser.add_argument(
        "-B",
        "--show-body",
        action="store_true",
        help="""
            Display the received body
        """,
    )

    args = parser.parse_args()
    run(
        address=args.bind_address,
        port=args.port,
        show_body=args.show_body,
    )
