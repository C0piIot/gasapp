from django.core.management.base import BaseCommand
from django.contrib.gis.geos import Point
from stations.models import Station
import urllib.request, json
from django.db import transaction

class Command(BaseCommand):
    help = 'Update stations pricing'
    json_url = 'https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/'
    
    def handle(self, *args, **options):
        with urllib.request.urlopen(self.json_url) as response:
            stations = json.loads(response.read())['ListaEESSPrecio']
            keys = {}
            with transaction.atomic():
                l = 0
                com = 0
                biodie = 0
                etan = 0
                gasonu =0
                gases =0
                prem95 = 0
                prem98 = 0
                for station in stations:
                    petrol95 = station['Precio Gasolina 95 E5'] or station['Precio Gasolina 95 E5 Premium']
                    petrol98 = station['Precio Gasolina 98 E5'] or station['Precio Gasolina 98 E10']
                    gasoil = station['Precio Gasoleo A'] or station['Precio Gasoleo Premium']
                    glp = station['Precio Gases licuados del petróleo']

                    if petrol98 or petrol95 or gasoil:

                        Station.objects.update_or_create(pk=station['IDEESS'],
                            defaults={
                                'pk': station['IDEESS'],
                                'name': station['Rótulo'].title(),
                                'postal_code': station['C.P.'],
                                'address': station['Dirección'].title(),
                                'opening_hours': station['Horario'],
                                'town': station['Localidad'].title(),
                                'city': station['Municipio'],
                                'state': station['Provincia'].title(),
                                'petrol95': petrol95.replace(',', '.') if petrol95 else None,
                                'petrol98': petrol98.replace(',', '.') if petrol98 else None,
                                'gasoil': gasoil.replace(',', '.') if gasoil else None,
                                'glp': glp.replace(',', '.') if glp else None,
                                'location': Point(
                                    float(station['Longitud (WGS84)'].replace(',','.')), 
                                    float(station['Latitud'].replace(',','.'))
                                )
                        })
