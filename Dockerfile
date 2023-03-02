FROM python:3-slim AS base
ENV PYTHONUNBUFFERED 1
ENV PYTHONDONTWRITEBYTECODE 1
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y python3-gdal libsqlite3-mod-spatialite python3-dev build-essential sqlite3 && \
    apt-get autoremove --purge -y
RUN pip install --upgrade pip
RUN mkdir /app
RUN mkdir /cache
WORKDIR /app
COPY requirements.txt /app/
RUN	pip install -r requirements.txt
CMD ["python", "manage.py", "runserver", "0.0.0.0:80"]

FROM base AS prod
COPY . /app/
RUN python manage.py collectstatic --no-input
CMD ["bash", "/app/entrypoint.sh"]

