FROM golang:1.14.2-buster AS gobuild

# run dependencies
RUN apt-get update \
    && apt-get install -y --no-install-recommends g++ make gcc git build-essential ca-certificates && \
    update-ca-certificates

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
RUN go build -a --ldflags '-extldflags "-static"' entrypoints/main.go


# copy executable file and certs to a pure container
FROM debian:buster

WORKDIR /app

COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
COPY --from=gobuild /goapp/main /app/go-graphql-srv

RUN chmod +rx -R /app && \
    adduser -S laisky
USER laisky

ENTRYPOINT [ "./go-graphql-srv" ]
CMD [ "--debug", "--addr=127.0.0.1:8080", "--dbaddr=127.0.0.1:27017" ]
