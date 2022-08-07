# Install

Gremlins can be installed via pre-compiled binaries or from source.

## Pre compiled binaries

### :material-linux: Linux

We don't have public repositories yet. To install, you have to download the package appropriate to your architecture/OS
and install it "manually".

=== ":material-debian: deb"

    Download a `.deb` file appropriate for your ARCH from
    the [release page](https://github.com/go-gremlins/gremlins/releases/latest), then install with:

    ```sh
    dpkg -i gremlins_{{ release.full_version }}_linux_amd64.deb
    ```

=== ":material-redhat: rpm"

    Download a `.rpm` file appropriate for your ARCH from
    the [release page](https://github.com/go-gremlins/gremlins/releases/latest), then install with:

    ```sh
    rpm -i gremlins_{{ release.full_version }}_linux_amd64.rpm
    ```

### :material-apple: MacOS

On macOS, you can use [Homebrew](https://brew.sh/). As of now, Gremlins uses only an Homebrew _tap_.

To install, you have to first _tap_ Gremlins' repository:

```sh
brew tap go-gremlins/tap #(1)
```

1. Doing this, your tap will refer directly to the Gremlins' _tap formula_ on GitHub. You can delete the _tap_ by
   "untapping" it:
   ```sh
   brew untap go-gremlins/tap
   ```

Then you can install it:

```sh
brew install gremlins
```

### :material-microsoft-windows: Windows

As of now, only manual installation is supported.
Download the appropriate release package from
the [release page](https://github.com/go-gremlins/gremlins/releases/latest),
extract the zip archive and copy the `.exe` file somewhere in your execution `PATH`.

### :material-docker: Docker

You can also run Gremlins using the official Docker image:

```shell
docker run --rm -v $(pwd):/app -w /app gogremlins/gremlins gremlins unleash .
```

### :material-bash: Manual install

Alternatively, you can download the binary for your OS/ARCH, _untar_ it.

For example, on GNU/Linux it could be:

```shell
tar -xvf gremlins_{{ release.full_version }}_linux_amd64.tar.gz
```

then copy it somewhere in `PATH`:

```shell
sudo cp gremlins_{{ release.full_version }}_linux_amd64/gremlins /usr/bin
```

## From source

### :fontawesome-brands-golang: Go install

Gremlins can be installed with the Go install command. Only the [Go compiler](https://go.dev) is needed.

```sh
go install https://github.com/go-gremlins/gremlins/cmd/gremlins@v{{ release.full_version }}
```

### :material-ninja: Ninja style

To build Gremlins you need the [Go compiler](https://go.dev), `make` and [golangci-lint](https://golangci-lint.run) for
linting. You can clone download the source tarball from
the [release page](https://github.com/go-gremlins/gremlins/releases/latest), then:

```sh
tar -xvf gremlins-{{ release.full_version }}.tar.gz
```

Ad then:

```sh
cd gremlins-{{ release.full_version }}
```

```sh
make
```

At this point, you can move the generated binary executable to a location of your choice.
