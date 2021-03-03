from django.views.generic.list import BaseListView
from django.contrib.gis.geos import Polygon, Point
from django.contrib.gis.db.models.functions import Distance
from django.http import JsonResponse
from .models import *

class StationsView(BaseListView):

    model = Station
    paginate_by = 100
    
    def get_queryset(self):
        queryset = super().get_queryset()
        if 'center' in self.request.GET:
            lat, lng = self.request.GET.get('center').split(',')
            p = Point(float(lat), float(lng), srid=4326)
            queryset = queryset.annotate(distance=Distance("location", p))
            queryset = queryset.filter(location__dwithin=(p, 0.4)).order_by('distance')
        return queryset.values(
            'pk', 'name', 'petrol95', 'petrol98', 'gasoil', 'address', 'city', 'postal_code', 'location', 'updated'
        )

    def render_to_response(self, context):
        return JsonResponse({ 'stations': [[
                s['pk'],
                s['name'],
                float(s['petrol95']) if s['petrol95'] else None,
                float(s['petrol98']) if s['petrol98'] else None,
                float(s['gasoil']) if s['gasoil'] else None,
                s['address'],
                s['city'],
                s['postal_code'],
                [coordinate for coordinate in s['location']],
                s['updated'].timestamp()
            ] for s in context['object_list']]})
