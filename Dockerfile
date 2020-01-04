FROM golang:1.13
WORKDIR /go/src/github.com/KageShiron/aspandoc
ENV GO111MODULE=on
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM kageshiron/pandoc
WORKDIR /root/
COPY --from=0 /go/src/github.com/KageShiron/aspandoc/app .
CMD ["./app"]
EXPOSE 8080
