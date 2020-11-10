# skynet-portal-rename

This script renames files on a [Skynet
Webportal](https://github.com/NebulousLabs/skynet-webportal) and then deletes
all the old empty directories. It will write the new directory names to disk for
reference as well as ensure that there are `.siadir` files in all the newly
created directories.

## Usage

There are two constants, `dirDepth` and `dirLength` that can be modified to
generate different desired file structures.

This script can also be used to clean up old attempts at renaming. It has
a `validDirStructure` function that will skip a rename if the file already is in
a directory that satisfies the expected filesystem structure defined by
`dirDepth` and `dirLength`. This also means that you can run this script
multiple times with incurring rework.

Lastly, you can take advantage of the empty directory deletion in the script by
supplying `delete-only` as an argument. This will skip the renaming and just
iterate over the filesystem and delete old empty directories.

