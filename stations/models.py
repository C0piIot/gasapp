from django.db import models
from django.contrib.gis.db import models as gis_models
from django.utils.translation import gettext_lazy as _

class Station(models.Model):
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
    location = gis_models.PointField(_('location'))

    class Meta:
        verbose_name = _('station')
        verbose_name_plural = _('stations')



    '''
https://gis.stackexchange.com/questions/141533/geodjango-find-all-points-within-radius
{'C.P.': '01240', 
'Dirección': 'CL MANISITU, 9',
 'Horario': 'L-D: 24H',

 'Latitud': '42,846028', 
 'Localidad': 'ALEGRIA-DULANTZI', 
 'Longitud (WGS84)': '-2,509361', 
 'Margen': 'D', 
 'Municipio': 'Alegría-Dulantzi',
  'Precio Biodiesel': None, 
  'Precio Bioetanol': None, 
  'Precio Gas Natural Comprimido': None, 
  'Precio Gas Natural Licuado': None,
   'Precio Gases licuados del petróleo': None, 
   'Precio Gasoleo A': '1,269',
    'Precio Gasoleo B': '0,726', 
    'Precio Gasolina 95 Protección': None,
     'Precio Gasolina  98': None, 
     'Precio Nuevo Gasoleo A': None, 
 'Provincia': 'ÁLAVA',
  'Remisión': 'dm', 
  'Rótulo': 'PREMIRA ENERGIA NORTE, S.L.', 
  'Tipo Venta': 'P', '% BioEtanol': '0,0', '% Éster metílico': '0,0',
   'IDEESS': '9381', 'IDMunicipio': '1', 'IDProvincia': '01', 'IDCCAA': '16'}'''

