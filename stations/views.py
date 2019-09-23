from django.views.generic.list import BaseListView
from django.contrib.gis.geos import Polygon, Point
from django.contrib.gis.db.models.functions import Distance
from django.http import JsonResponse
from .models import *

class StationsView(BaseListView):

    model = Station
    paginate_by = 300

    def get_queryset(self):
        queryset = super().get_queryset()
        if 'center' in self.request.GET:
            lat, lng = self.request.GET.get('center').split(',')
            queryset = queryset.annotate(distance=Distance("location", Point(float(lat), float(lng), srid=4326))).order_by('distance')
        return queryset

    def render_to_response(self, context):
        return JsonResponse({ 'stations': [[
                s.pk,
                s.name,
                s.petrol95,
                s.petrol98,
                s.gasoil,
                s.address,
                s.town,
                s.city,
                s.postal_code,
                [coordinate for coordinate in s.location]
            ] for s in context['object_list']]})