FROM golang:1.8-alpine

RUN apk --no-cache add -t build-deps build-base go git \
	&& apk --no-cache add ca-certificates

WORKDIR /go/src/app
COPY ./src/ .

RUN go-wrapper download
RUN go-wrapper install

RUN mkdir -p ~/.config/itomych/calendar-bot && mkdir ~/.credentials
COPY config.yaml /root/.config/itomych/calendar-bot/
COPY itomych-calendar-bot.json /root/.credentials/

CMD ["go-wrapper", "run"]
