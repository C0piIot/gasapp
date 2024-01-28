FROM python:3.11-slim AS base
ENV PYTHONUNBUFFERED 1
ENV PYTHONDONTWRITEBYTECODE 1
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y --no-install-recommends python3-gdal libsqlite3-mod-spatialite python3-dev build-essential sqlite3 yarnpkg && \
    apt-get autoremove --purge -y
RUN pip install --upgrade pip
RUN mkdir /app
RUN mkdir /cache
WORKDIR /app
COPY requirements.txt package.json yarn.lock /app/
RUN	pip install -r requirements.txt
RUN yarnpkg install
CMD ["python", "manage.py", "runserver", "0.0.0.0:80"]

FROM base AS prod
ARG DEBUG False
ARG SENTRY_DSN=https://public@sentry.example.com/1
COPY . /app/
RUN python manage.py collectstatic --no-input
RUN python manage.py compress
CMD ["bash", "/app/entrypoint.sh"]

