FROM golang:1.25-alpine AS apiBuilder
WORKDIR /opt
RUN apk update && apk add --no-cache make
COPY . /opt/
RUN go mod download
ARG GIT_TAG=?
RUN make build GIT_TAG=${GIT_TAG}

FROM alpine
ARG ENVIRONMENT=production
WORKDIR /opt
RUN apk update && apk add --no-cache curl ca-certificates ffmpeg-dev
COPY --from=apiBuilder /opt/k0pern1cus /opt/k0pern1cus
EXPOSE 8080
RUN ulimit -n 100000
CMD [ "./k0pern1cus" ]
