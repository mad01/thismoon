FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY tools/suspenders ./tools/suspenders
RUN CGO_ENABLED=0 GOOS=linux go build -o /suspenders ./tools/suspenders/cmd/suspenders/

FROM alpine:3.21
RUN apk add --no-cache bash git
RUN addgroup -S testgroup && adduser -S testuser -G testgroup
COPY --from=builder /suspenders /usr/local/bin/suspenders
USER testuser
WORKDIR /home/testuser
RUN git config --global user.email "test@test.com" && \
    git config --global user.name "Test User"
ENTRYPOINT ["/usr/local/bin/suspenders"]
