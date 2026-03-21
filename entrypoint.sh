#!/bin/bash

python manage.py migrate
sqlite3 /app/db.sqlite3 < /app/pragma.sql

while true; do python manage.py update_prices; echo "Prices updated"; sleep 3600; done&
uwsgi /app/uwsgi.ini
