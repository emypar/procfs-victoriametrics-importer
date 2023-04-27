#! /usr/bin/env python3

# Capture a snapshot of /proc filesystem

import argparse
import json
import os
import pwd
import re
import shutil
import sys
import time
from typing import List, Optional, Set, Tuple, Union

DEFAULT_PROC_DIR = "/proc"
uid = os.getuid()
DEFAULT_DST_DIR = os.path.join(
    (f"/tmp/{pwd.getpwuid(uid).pw_name}" if uid != 0 else "/var/proc-snap"),
    "snap",
    "%FT%T%z",
    "proc",
)
DEFAULT_REPEAT = None
DEFAULT_PAUSE_SEC = 10
SingleMatch = Union[str, re.Pattern, Set[str]]
Match = Union[SingleMatch, List[SingleMatch]]
IgnoreList = List[Tuple[Match, Match]]

METADATA_FILE = ".metadata.json"

# All paths below are relative to proc root:
IGNORE_PATH_LIST = [
    # /proc/PID and /proc/PID/task/TID exceptions:
    (
        re.compile("\d+(/task/\d+)?"),
        set(
            [
                "attr",
                "clear_refs",
                "map_files",
                "mem",
                # "net",
                "ns",
                "pagemap",
                "smaps_rollup",
                "stack",
            ]
        ),
    ),
    # /proc:
    (
        "",
        set(
            [
                "bus",
                "clear_refs",
                "config.gz",
                "driver",
                "ide",
                "kallsyms",
                "kcore",
                "keys",
                "kmsg",
                "ksyms",
                "malloc",
                "modules",
                "mttr",
                "pagemap",
                "pci",
                "scsi",
                "smaps_rollup",
                "sys",
                "sysrq-trigger",
                "sysvipc",
                "tty",
            ]
        ),
    ),
]


def single_match(match: SingleMatch, val: str) -> bool:
    if isinstance(match, str):
        return val == match
    elif isinstance(match, re.Pattern):
        return match.match(val) is not None
    elif isinstance(match, set):
        return val in match
    else:
        return False


def is_ignored(dirpath: str, name: str) -> bool:
    return any(
        single_match(m[0], dirpath) and single_match(m[1], name)
        for m in IGNORE_PATH_LIST
    )


def make_ignore(root: str = DEFAULT_PROC_DIR):
    root = os.path.normpath(root)
    root_slash = root + "/"
    skip_root_index = len(root_slash)

    def ignore(dirpath: str, names: List[str]) -> Set[str]:
        if dirpath == root:
            dirpath = ""
        elif dirpath.startswith(root_slash):
            dirpath = dirpath[skip_root_index:]
        return set(name for name in names if is_ignored(dirpath, name))

    return ignore


def do_snap(proc_root, dst_root):
    start_time = time.time()

    # Use copy instead of copy2 to get the timestamp when the snap was created.
    try:
        shutil.copytree(
            proc_root,
            dst_root,
            copy_function=shutil.copy,
            symlinks=True,
            ignore=make_ignore(proc_root),
            ignore_dangling_symlinks=True,
            dirs_exist_ok=True,
        )
    except (shutil.Error) as e:
        for _, _, errormsg in e.args[0]:
            print(errormsg, file=sys.stderr)
    d_time = time.time() - start_time
    print(
        f"{proc_root!r} snapped to {dst_root!r} in {d_time:.02f} sec", file=sys.stderr
    )


def do_post_snap_fixes(dst_root, user: Optional[str] = None):
    self_path = os.path.join(dst_root, "self")
    thread_self_path = os.path.join(dst_root, "thread-self")
    try:
        os.unlink(self_path)
        os.symlink("1", self_path)
        os.unlink(thread_self_path)
        os.symlink("1/task/1", thread_self_path)
        os.system(f"chmod -R a+r-w {dst_root}")
        if user is not None:
            os.system(f"chown -R {user} {dst_root}")
    except KeyboardInterrupt as e:
        raise e
    except Exception as e:
        print(e, file=sys.stderr)


def add_metadata(dst_root):
    uname_info = os.uname()
    clktck = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
    with open(os.path.join(dst_root, METADATA_FILE), "wt") as f:
        json.dump(
            {
                "arch": uname_info.machine,
                "os": uname_info.sysname,
                "release": uname_info.release,
                "version": uname_info.version,
                "clktck": clktck,
            },
            f,
            indent=2,
        )
        f.write("\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-p",
        "--proc-root",
        default=DEFAULT_PROC_DIR,
        help="""
            procfs root dir, default: %(default)s
        """,
    )
    parser.add_argument(
        "-d",
        "--destination",
        default=DEFAULT_DST_DIR,
        help="""
            Destination root dir passed through strftime, default: %(default)s
        """,
    )
    parser.add_argument(
        "-u",
        "--user",
        help="""
            Change snap ownership to USER.
        """,
    )

    parser.add_argument(
        "-r",
        "--repeat",
        type=int,
        default=DEFAULT_REPEAT,
        help="""
            Repeat the snap, default: %(default)s
        """,
    )
    parser.add_argument(
        "-s",
        "--pause",
        type=float,
        default=DEFAULT_PAUSE_SEC,
        help="""
            Pause between repeats in seconds, default: %(default)f
        """,
    )

    args = parser.parse_args()
    repeat = args.repeat or 1
    pause_sec = args.pause
    next_snap_ts = None
    for _ in range(repeat):
        if next_snap_ts is not None:
            pause_till_next_snap = next_snap_ts - time.time()
            if pause_till_next_snap > 0:
                time.sleep(pause_till_next_snap)
                next_snap_ts += pause_sec
        else:
            next_snap_ts = time.time() + pause_sec
        dst_root = time.strftime(args.destination)
        do_snap(args.proc_root, dst_root)
        do_post_snap_fixes(dst_root, user=args.user)
        add_metadata(dst_root)
