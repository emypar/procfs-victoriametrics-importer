#! /bin/bash

# Apply job code formatting tools:

case "$0" in
    /*|*/*) this_dir=$(cd $(dirname $0) && pwd);;
    *) this_dir=$(cd $(dirname $(which $0)) && pwd);;
esac

set -e
cd $this_dir
set -x
autoflake --in-place --remove-all-unused-imports --ignore-init-module-imports --recursive .
isort .
black .

