# Install

go-rae-gremlins publishes pre-compiled binaries as assets of its GitHub releases. You can also install it from source.

## Pre-compiled binaries

Download the archive for your OS and architecture from the
[release page](https://github.com/OpenRAE/go-rae-gremlins/releases/latest). Each release has archives for Linux
(386, amd64, arm64), macOS (amd64, arm64), and Windows (386, amd64, arm64), and a `checksums.txt` file.

Extract the archive and copy the `gremlins` binary to a directory in your `PATH`. For example, on GNU/Linux:

```shell
tar -xvf gremlins_<version>_linux_amd64.tar.gz
sudo cp gremlins /usr/local/bin
```

## From source

### Go install

Only the [Go compiler](https://go.dev) is necessary:

```sh
go install github.com/OpenRAE/go-rae-gremlins/cmd/gremlins@v<version>
```

### Build

To build Gremlins, you need the [Go compiler](https://go.dev) and `make`:

```sh
git clone https://github.com/OpenRAE/go-rae-gremlins.git
cd go-rae-gremlins
make build
```

The binary is `dist/bin/gremlins`.
