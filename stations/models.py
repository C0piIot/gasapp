from django.db import models, transaction
from django.contrib.gis.db import models as gis_models
from django.utils.translation import gettext_lazy as _
from django.contrib.gis.geos import Point
from urllib import request
from xml.etree import cElementTree as ET

ns                  = '{http://schemas.datacontract.org/2004/07/ServiciosCarburantes}%s'
STATION             = ns % 'EESSPrecio'
PETROL95            = ns % 'Precio_x0020_Gasolina_x0020_95_x0020_E5'
PETROL95_ALTERNATE  = ns % 'Precio_x0020_Gasolina_x0020_95_x0020_E5_x0020_Premium'
PETROL98            = ns % 'Precio_x0020_Gasolina_x0020_98_x0020_E5'
PETROL98_ALTERNATE  = ns % 'Precio_x0020_Gasolina_x0020_98_x0020_E10'
GASOIL              = ns % 'Precio_x0020_Gasoleo_x0020_A'
GASOIL_ALTERNATE    = ns % 'Precio_x0020_Gasoleo_x0020_Premium'
GLP                 = ns % 'Precio_x0020_Gases_x0020_licuados_x0020_del_x0020_petróleo'
ID                  = ns % 'IDEESS'
NAME                = ns % 'Rótulo'
POSTAL_CODE         = ns %'C.P.'
ADDRESS             = ns % 'Dirección'
OPENING_HOURS       = ns % 'Horario'
TOWN                = ns %'Localidad'
CITY                = ns % 'Municipio'
STATE               = ns %'Provincia'
LONG                = ns % 'Longitud_x0020__x0028_WGS84_x0029_'
LAT                 = ns % 'Latitud'


class Station(models.Model):
    PRICES_URL = 'https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/'

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
        with request.urlopen(request.Request(cls.PRICES_URL, headers={'Accept': 'application/xml'})) as response:
            with transaction.atomic():
                for event, elem in ET.iterparse(response, events=("end",)):
                    if event == 'end' and elem.tag == STATION:
                        cls.update_station(elem)
    
    @classmethod
    def update_station(cls, elem):
        petrol95 = elem.find(PETROL95).text or elem.find(PETROL95_ALTERNATE).text
        petrol98 = elem.find(PETROL98).text or elem.find(PETROL98_ALTERNATE).text
        gasoil = elem.find(GASOIL).text or elem.find(GASOIL_ALTERNATE).text
        glp = elem.find(GLP).text

        if petrol95 or petrol98 or gasoil or glp:
            cls.objects.update_or_create(pk=elem.find(ID).text,
                defaults={
                    'pk': elem.find(ID).text,
                    'name': elem.find(NAME).text.title(),
                    'postal_code': elem.find(POSTAL_CODE).text,
                    'address': elem.find(ADDRESS).text.title(),
                    'opening_hours': elem.find(OPENING_HOURS).text,
                    'town': elem.find(TOWN).text.title(),
                    'city': elem.find(CITY).text,
                    'state': elem.find(STATE).text.title(),
                    'petrol95': petrol95.replace(',', '.') if petrol95 else None,
                    'petrol98': petrol98.replace(',', '.') if petrol98 else None,
                    'gasoil': gasoil.replace(',', '.') if gasoil else None,
                    'glp': glp.replace(',', '.') if glp else None,
                    'location': Point(
                        float(elem.find(LONG).text.replace(',','.')), 
                        float(elem.find(LAT).text.replace(',','.'))
                    )
            })
