#!/bin/bash

python manage.py migrate
while true; do python manage.py update_prices; echo "Prices updated"; sleep 3600; done&
uwsgi /code/docker/uwsgi.ini
