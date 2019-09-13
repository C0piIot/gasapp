FROM python:alpine
ENV PYTHONUNBUFFERED 1
RUN mkdir /code
WORKDIR /code
COPY requirements.txt /code/
RUN apk update && \
    apk --no-cache upgrade && \
    apk add --no-cache \
    	--repository http://dl-cdn.alpinelinux.org/alpine/edge/testing \
  		--repository http://dl-cdn.alpinelinux.org/alpine/edge/main \
  		gdal geos sqlite libspatialite libspatialite-dev proj && \
  	apk add \
  		--repository http://dl-cdn.alpinelinux.org/alpine/edge/testing \
  		--repository http://dl-cdn.alpinelinux.org/alpine/edge/main \
  		--no-cache --virtual .build-deps gcc python3-dev gdal-dev build-base && \
	pip install --upgrade pip && \
	pip install -r requirements.txt && \
	apk del --no-cache .build-deps && \
  ln -s /usr/lib/libproj.so.15 /usr/lib/libproj.so

#apk add --no-cache --virtual .build-deps postgresql-dev gcc python3-dev musl-dev linux-headers 