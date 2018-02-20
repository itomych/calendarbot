# CalendarBot
>A simple bot that manages shared Google Calendar

We need a bot that would manage our GCal user for a meeting room. The bot checks GCal with some configurable interval and 
accepts or declines upcoming events depending on the conflicts

# Installation

1. Follow the [Google Calendar API Go guide](https://developers.google.com/google-apps/calendar/quickstart/go) to setup your project and get credetials.
   Save the credentials to `client_credentials.json` in `src` folder.

1. Create `config.yaml` in the root folder like this sample:
```yaml
email: meetingroom@mydomain.com
checkinterval: 60
```
1. Build Docker container `docker build -t itomychstudio/calendarbot .`

1. First run should be done in the interactive mode to get the token from Google OAuth2 `docker run -it --name calendarbot itomychstudio/calendarbot:latest`

1. Afterwards, you can run the container in the detached mode `docker run -d --name calendarbot --restart always itomychstudio/calendarbot:latest`.

If you cannot run the container in the interactive mode, run the app locally and copy the ~/.credentials/itomych-calendar-bot.json file to the root folder of the project. 
It will be automatically copied to the container on docker build.