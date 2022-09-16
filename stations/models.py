from django.db import models, transaction
from django.contrib.gis.db import models as gis_models
from django.utils.translation import gettext_lazy as _
from django.contrib.gis.geos import Point
import urllib.request, json


class Station(models.Model):
    json_url = 'https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/'

    name = models.CharField(max_length=200)
    updated = models.DateTimeField(auto_now=True)
    postal_code = models.CharField(_('postal code'), max_length=8)
    address = models.CharField(_('address'), max_length=200)
    opening_hours = models.CharField(_('opening hours'), max_length=200)
    town = models.CharField(_('town'), max_length=200)
    city = models.CharField(_('city'), max_length=200)
    state = models.CharField(_('state'), max_length=200)
    gasoil = models.DecimalField(_('gasoil'), max_digits=6, decimal_places=3, blank=True, null=True)
    petrol95 = models.DecimalField(_('gasolina 95'), max_digits=6, decimal_places=3, blank=True, null=True)
    petrol98 = models.DecimalField(_('gasolina 98'), max_digits=6, decimal_places=3, blank=True, null=True)
    glp = models.DecimalField(_('GLP'), max_digits=6, decimal_places=3, blank=True, null=True)
    location = gis_models.PointField(_('location'))

    class Meta:
        verbose_name = _('station')
        verbose_name_plural = _('stations')

    
    @classmethod
    def update_prices(cls):
        with urllib.request.urlopen(cls.json_url) as response:
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

                        cls.objects.update_or_create(pk=station['IDEESS'],
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

