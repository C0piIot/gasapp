FROM python:alpine
ENV PYTHONUNBUFFERED 1
RUN mkdir /code
WORKDIR /code
COPY requirements.txt /code/
RUN apk update && \
    apk --no-cache upgrade && \
    apk add gdal && \
    pip install --upgrade pip && \
    pip install -r requirements.txt