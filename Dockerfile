FROM pypy:3-slim AS base
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y --no-install-recommends gdal-bin libgdal-dev libgeos-c1v5 libsqlite3-mod-spatialite build-essential sqlite3 yarnpkg && \
    apt-get autoremove --purge -y
RUN pip install --upgrade pip
RUN mkdir /app
RUN mkdir /cache
WORKDIR /app
COPY requirements.txt package.json yarn.lock /app/
RUN	pip install -r requirements.txt
RUN yarnpkg install
CMD ["pypy3", "manage.py", "runserver", "0.0.0.0:80"]
ARG BUILD_VERSION=dev
ENV BUILD_VERSION=$BUILD_VERSION
ARG GIT_REV=HEAD
ENV GIT_REV=$GIT_REV

FROM base AS prod
ARG DEBUG False
COPY . /app/
RUN pypy3 manage.py collectstatic --no-input
RUN pypy3 manage.py compress
CMD ["bash", "/app/entrypoint.sh"]

