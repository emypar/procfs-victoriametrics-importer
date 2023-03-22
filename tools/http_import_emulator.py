#! /usr/bin/env python3

# Simulate HTTP import endpoint: accept and display requests.

import argparse
import gzip
import http.client
import http.server
import re
import socketserver
from http import HTTPStatus
from typing import List, Optional, Tuple, Union

DEFAULT_PORT = 8080

ChunkExtension = Union[str, Tuple[str, str]]


def parse_chunk_size_and_extension(
    chunk_sz_ext: str,
) -> Tuple[int, List[ChunkExtension]]:
    sz_ext = re.split(r"\s*;\s*", chunk_sz_ext)
    return int(sz_ext[0], base=16), [
        re.split(r"\s*=\s*", e.strip()) for e in sz_ext[1:]
    ]


class HTTPRequestHandler(http.server.BaseHTTPRequestHandler):
    def read_chunk(self) -> Optional[bytes]:
        chunk_sz_ext = b""
        # Read until '\r':
        while True:
            b = self.rfile.read(1)
            if len(b) == 0:
                return None
            if b == b"\r":
                break
            chunk_sz_ext += b
        # Expect '\n':
        b = self.rfile.read(1)
        if b != b"\n":
            return None
        # Parse chunk_sz_ext:
        chunk_sz, chunk_ext = parse_chunk_size_and_extension(str(chunk_sz_ext, "utf-8"))
        self.log_message(
            "chunk size=%d, chunk extensions=%s", chunk_sz, repr(chunk_ext)
        )
        # Read the actual chunk:
        if chunk_sz > 0:
            chunk = self.rfile.read(chunk_sz)
        else:
            chunk = b""
        if len(chunk) != chunk_sz:
            return None
        # Verify '\r\n' after the chunk:
        if self.rfile.read(2) != b"\r\n":
            return None
        return chunk

    def do_POST(self):
        self.log_message("%s\n%s", self.requestline, self.headers)

        content_encoding = self.headers.get("Content-Encoding")
        if content_encoding not in {None, "gzip"}:
            self.send_error(
                HTTPStatus.UNSUPPORTED_MEDIA_TYPE,
                HTTPStatus.UNSUPPORTED_MEDIA_TYPE.description,
                explain=f"Content-Encoding: {content_encoding} not supported"
            )
            return

        content_type = (
            self.headers.get_content_type() or self.headers.get_default_type()
        )
        content_main_type, content_sub_type = content_type.split("/")
        if content_main_type != "text":
            self.send_error(
                HTTPStatus.NOT_IMPLEMENTED, 
                message=HTTPStatus.NOT_IMPLEMENTED.description,
                explain=f"Content-Type: {content_type} not supported"
            )
            return

        transfer_encoding = self.headers.get("Transfer-Encoding")
        if transfer_encoding == "chunked":
            body = b""
            if self.headers.get("Expect") == "100-continue":
                self.send_response(HTTPStatus.CONTINUE)
                self.end_headers()
            while True:
                chunk = self.read_chunk()
                if chunk is None:
                    self.send_error(
                        HTTPStatus.UNPROCESSABLE_ENTITY,
                        HTTPStatus.UNPROCESSABLE_ENTITY.description,
                        explain="Invalid chunk"
                    )
                    return
                if len(chunk) == 0:
                    break
                body += chunk
        elif transfer_encoding is None:
            try:
                content_length = int(self.headers["Content-Length"])
            except (ValueError, TypeError, KeyError):
                self.send_error(
                    HTTPStatus.LENGTH_REQUIRED, 
                    HTTPStatus.LENGTH_REQUIRED.description,
                    explain="missing Content-Length"
                )
                return
            body = self.rfile.read(content_length)
            if len(body) != content_length:
                self.send_error(
                    HTTPStatus.UNPROCESSABLE_ENTITY,
                    HTTPStatus.UNPROCESSABLE_ENTITY.description,
                )
                return
        else:
            self.send_error(
                HTTPStatus.UNPROCESSABLE_ENTITY,
                message=HTTPStatus.UNPROCESSABLE_ENTITY.description,
                explain=f"Transfer-Encoding: {transfer_encoding} not supported"
            )

        if content_encoding == "gzip":
            body = gzip.decompress(body)
        self.log_message("body:\n%s", str(body, "utf-8"))
        self.send_response(HTTPStatus.OK)
        self.flush_headers()

    def do_GET(self):
        self.log_message(f""""{self.requestline}"\n{self.headers}""")
        self.send_response(HTTPStatus.OK)

    do_HEAD = do_GET
    do_PUT = do_POST


def run(
    address="",
    port=DEFAULT_PORT,
):
    with socketserver.ThreadingTCPServer((address, port), HTTPRequestHandler) as httpd:
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
    args = parser.parse_args()
    run(address=args.bind_address, port=args.port)
