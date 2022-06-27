all: app

app: *.go go.mod go.sum
	GOOS=linux GOARCH=amd64 go build -o app
