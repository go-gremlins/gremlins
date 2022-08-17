# Docker

Gremlins can be used in CI pipelines using the prebuilt Docker images.

## As a pipeline step

In the _continuous integration_ tool of your choice, you can execute a step using the following syntax:

```shell
docker run --rm -v $(pwd):/app -w /app gogremlins/gremlins:{{ release.full_version }} gremlins unleash .
```

The exact way to specify a runner step in the pipeline script depends on the tool of choice.

## As a stage in the Dockerfile

Gremlins can be also run as a stage in the `Dockerfile`.

```dockerfile
FROM gogremlins/gremlins:{{ release.full_version }} AS mutation-testing
WORKDIR /my/project/source
RUN gremlins unleash
```

For further details, please refer to
the [Docker multi stage builds documentation](https://docs.docker.com/develop/develop-images/multistage-build/).