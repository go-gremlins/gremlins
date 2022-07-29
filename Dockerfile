FROM golang:1.18

COPY gremlins /usr/bin
RUN adduser --disabled-password --gecos "" nonroot
USER nonroot:nonroot

CMD ["gremlins"]