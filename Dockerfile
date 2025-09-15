FROM golang:1.25 AS apiBuilder
WORKDIR /opt
RUN apt-get update && apt-get install -y make
COPY . /opt/
RUN go mod download
ARG GIT_TAG=?
RUN make build GIT_TAG=${GIT_TAG}

FROM ubuntu:22.04
ARG ENVIRONMENT=production
WORKDIR /opt
RUN apt-get update && apt-get install -y curl ca-certificates ffmpeg && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
COPY --from=apiBuilder /opt/k0pern1cus /opt/k0pern1cus
EXPOSE 8080
RUN ulimit -n 100000
CMD [ "./k0pern1cus" ]