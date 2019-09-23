FROM python
ENV PYTHONUNBUFFERED 1
RUN mkdir /code
WORKDIR /code
COPY requirements.txt /code/
RUN apt-get -y update && \
    apt-get -y dist-upgrade && \
    apt-get -y install binutils libproj-dev gdal-bin && \
	   pip install --upgrade pip && \
	   pip install -r requirements.txt