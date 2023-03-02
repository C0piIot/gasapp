FROM python:3-slim
ENV PYTHONUNBUFFERED 1
ENV PYTHONDONTWRITEBYTECODE 1
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y python3-gdal libsqlite3-mod-spatialite python3-dev build-essential sqlite3 && \
    apt-get autoremove --purge -y
RUN pip install --upgrade pip
RUN mkdir /code
RUN mkdir /cache
WORKDIR /code
COPY requirements.txt /code/
RUN	pip install -r requirements.txt
COPY . /code/
RUN python manage.py collectstatic --no-input
CMD ["bash", "/code/docker/entrypoint.sh"]
#CMD ["python", "manage.py", "runserver", "0.0.0.0:80"]