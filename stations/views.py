from django.views.generic.list import BaseListView
from django.http import JsonResponse
from .models import *

class StationsView(BaseListView):

    model = Station
    paginate_by = 100

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