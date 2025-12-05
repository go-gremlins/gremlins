FROM golang:1.25

COPY gremlins /usr/bin
RUN adduser --disabled-password --gecos "" nonroot
USER nonroot:nonroot

CMD ["gremlins"]