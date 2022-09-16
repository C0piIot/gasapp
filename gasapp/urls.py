from django.urls import path
from django.views.generic.base import TemplateView
from stations.views import *
from django.views.decorators.cache import cache_page

urlpatterns = [
    path('', cache_page(60 * 60 * 24)(TemplateView.as_view(template_name='home.html')), name='home'),
    path('stations/', cache_page(60 * 60 * 24)(StationsView.as_view()), name="stations"),
    path('offline/', cache_page(60 * 60 * 24)(TemplateView.as_view(template_name='offline.html')), name='offline'),
    path('update_prices/', cache_page(60 * 60)(UpdatePricesView.as_view()), name='update_prices')
]
