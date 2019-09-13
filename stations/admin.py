from django.contrib.gis import admin 
from .models import *
# Register your models here.
@admin.register(Station)
class StationAdmin(admin.OSMGeoAdmin):
	ate_hierarchy = 'updated'
	search_fields = ('name', 'town', 'city', 'address', 'postal_code',)
	list_display = ('name', 'updated', 'town', 'city', 'petrol95', 'petrol98', 'gasoil',)