#! /usr/bin/env python3

# Simulate HTTP import endpoint: accept and display requests.

import argparse
import gzip
import http.client
import http.server
import re
import socketserver
from http import HTTPStatus
from typing import BinaryIO, Callable, Optional

DEFAULT_PORT = 8080


class BodyError(Exception):
    pass


class BodyReader:
    def __init__(self, fh: BinaryIO, content_length: int = 0):
        if content_length <= 0:
            raise BodyError(f"{content_length}: Invalid Content-Length")
        self._fh = fh
        self._content_length = content_length
        self._n_read = 0

    def read(self, n: Optional[int]) -> bytes:
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


class ChunkedBodyReader:
    def __init__(self, fh: BinaryIO, fmt_logger: Optional[Callable]):
        self._fh = fh
        self._fmt_logger = fmt_logger
        self._chunk_sz, self._chunk_ext = None, None
        self._last_chunk = False

    def _parse_chunk_size_and_extension(self, chunk_sz_ext: bytes):
        try:
            sz_ext = re.split(r"\s*;\s*", str(chunk_sz_ext, "utf-8"))
            self._chunk_sz = int(sz_ext[0], base=16)
            self._chunk_ext = [re.split(r"\s*=\s*", e.strip()) for e in sz_ext[1:]]
        except (ValueError, TypeError, IndexError, UnicodeDecodeError):
            raise BodyError("Cannot process chunk header")

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
        self._parse_chunk_size_and_extension(chunk_sz_ext)
        if self._fmt_logger is not None:
            self._fmt_logger(
                "Chunk size: %d, extensions: %s",
                self._chunk_sz,
                repr(self._chunk_ext),
            )
        self._chunk_read = 0

    def read(self, n: Optional[int] = None) -> bytes:
        b = b""
        n_read = 0
        while (n is None or n_read < n) and not self._last_chunk:
            if self._chunk_sz is None:
                self._locate_next_chunk()
                if self._chunk_sz is None:
                    raise BodyError("Cannot locate chunk start")
            self._last_chunk = self._chunk_sz == 0
            if not self._last_chunk:
                to_read = min(n - n_read, self._chunk_sz - self._chunk_read)
                chunk_b = self._fh.read(to_read)
                if len(chunk_b) != to_read:
                    raise BodyError("Cannot read full chunk")
                if len(b) > 0:
                    b += chunk_b
                else:
                    b = chunk_b
                n_read += to_read
                self._chunk_read += to_read
            if self._chunk_read == self._chunk_sz:
                # Verify '\r\n' after the chunk:
                if self._fh.read(2) != b"\r\n":
                    raise BodyError("Cannot locate chunk end")
                # Done w/ this chunk:
                self._chunk_sz, self._chunk_ext = None, None
        return b


class DecodeGzip:
    def __init__(self, fh: BinaryIO):
        self._fh = gzip.open(fh, "rb")

    def read(self, n: Optional[int] = None) -> bytes:
        return self._fh.read(n)


SUPPORTED_ENCODING = {
    "gzip": DecodeGzip,
    "x-gzip": DecodeGzip,
}


class HTTPRequestHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.log_message("%s\n%s", self.requestline, self.headers)

        transfer_encodings = set(
            re.split(r"\s*,\s*", self.headers.get("Transfer-Encoding", "").strip())
        )
        if "chunked" in transfer_encodings:
            body_reader = ChunkedBodyReader(self.rfile, self.log_message)
            transfer_encodings.discard("chunked")
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
        for encoding in transfer_encodings:
            if encoding == "":
                pass
            elif encoding in SUPPORTED_ENCODING:
                body_reader = SUPPORTED_ENCODING[encoding](body_reader)
                break
            else:
                self.send_error(
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE,
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE.description,
                    explain=f"{encoding}: unsupported Transfer-Encoding",
                )
                return

        content_encodings = re.split(
            r"\s*,\s*", self.headers.get("Content-Encoding", "").strip()
        )
        for encoding in content_encodings:
            if encoding == "":
                pass
            elif encoding in SUPPORTED_ENCODING:
                body_reader = SUPPORTED_ENCODING[encoding](body_reader)
                break
            else:
                self.send_error(
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE,
                    HTTPStatus.UNSUPPORTED_MEDIA_TYPE.description,
                    explain=f"{encoding}: unsupported Content-Encoding",
                )
                return

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
