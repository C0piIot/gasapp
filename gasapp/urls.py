from django.contrib import admin
from django.urls import path
from django.views.generic.base import TemplateView
from stations.views import *

urlpatterns = [
    path('', TemplateView.as_view(template_name='home.html'), name='home'),
    path('stations/', StationsView.as_view(), name="stations"),
    path('admin/', admin.site.urls),
]
