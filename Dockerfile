FROM golang:1.9-alpine as build

RUN go version

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

RUN apk add --no-cache --update git &&\
    go get -u gopkg.in/alecthomas/gometalinter.v2 && \
    ln -s /go/bin/gometalinter.v2 /go/bin/gometalinter && \
    gometalinter --install --force

COPY . /go/src/github.com/itomych/calendarbot
WORKDIR /go/src/github.com/itomych/calendarbot

RUN cd src && go-wrapper download && go-wrapper install && go test -v $(go list -e ./... | grep -v vendor)

RUN gometalinter --disable-all --deadline=300s --vendor --enable=vet --enable=vetshadow --enable=golint \
    --enable=staticcheck --enable=ineffassign --enable=goconst --enable=errcheck --enable=unconvert \
    --enable=deadcode  --enable=gosimple --exclude=test --exclude=mock ./...

RUN go build -o calendarbot -ldflags "-X main.revision=$(git rev-parse --abbrev-ref HEAD)-$(git describe --abbrev=7 --always --tags)-$(date +%Y%m%d-%H:%M:%S) -s -w" ./src

FROM alpine:3.7

LABEL key="maitainer" value="Mikhail Merkulov <mikhail.m@itomy.ch>"

RUN apk add --update --no-cache tzdata
RUN apk add ca-certificates && rm -rf /var/cache/apk/*

WORKDIR /srv/

COPY --from=build /go/src/github.com/itomych/calendarbot/calendarbot /srv/

COPY init.sh /init.sh
RUN chmod +x /init.sh

RUN mkdir -p ~/.config/itomych/calendar-bot && mkdir ~/.credentials
COPY config.yaml /root/.config/itomych/calendar-bot/
COPY itomych-calendar-bot.json /root/.credentials/
COPY ./src/client_credentials.json /srv/

CMD ["/srv/calendarbot"]
ENTRYPOINT ["/init.sh"]