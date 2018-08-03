FROM umputun/baseimage:buildgo-latest as build

WORKDIR /go/src/github.com/itomych/calendarbot

ADD src /go/src/github.com/itomych/calendarbot/app
ADD .git /go/src/github.com/itomych/calendarbot/.git

RUN cd app && go test ./...

RUN gometalinter --disable-all --deadline=300s --vendor --enable=vet --enable=vetshadow --enable=golint \
    --enable=staticcheck --enable=ineffassign --enable=goconst --enable=errcheck --enable=unconvert \
    --enable=deadcode  --enable=gosimple --enable=gas --exclude=test --exclude=mock --exclude=vendor ./...

#RUN mkdir -p target && /script/coverage.sh

RUN \
    version=$(/script/git-rev.sh) && \
    echo "version $version" && \  
    go build -o calendarbot -ldflags "-X main.revision=${version} -s -w" ./app

FROM umputun/baseimage:app-latest

LABEL key="maitainer" value="Mikhail Merkulov <mikhail.m@itomy.ch>"

RUN apk add --update --no-cache tzdata
RUN apk add ca-certificates && rm -rf /var/cache/apk/*

WORKDIR /srv/

COPY --from=build /go/src/github.com/itomych/calendarbot/calendarbot /srv/

COPY init.sh /srv/init.sh
RUN chmod +x /srv/*.sh

RUN mkdir -p ~/.config/itomych/calendar-bot && mkdir ~/.credentials
COPY *.json /root/.credentials/
COPY ./src/client_credentials.json /srv/

CMD ["/srv/calendarbot"]
ENTRYPOINT ["/init.sh"]