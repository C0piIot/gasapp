from django.core.management.base import BaseCommand
import urllib.request, json

class Command(BaseCommand):
    help = 'Update stations pricing'
    json_url = 'https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/'
    
    def handle(self, *args, **options):
        with urllib.request.urlopen(self.json_url) as response:
            stations = json.loads(response.read())['ListaEESSPrecio']
            keys = {}
            for station in stations:
                for k in station:
                    if k in keys:
                        keys[k] += 1 if station[k] else 0
                    else:
                        keys[k] = 1 if station[k] else 0
            print(keys)

'''
'C.P.': 10341, 
'Dirección': 10341, 
'Horario': 10341, 
'Latitud': 10341, 
'Localidad': 10341, 
'Longitud (WGS84)': 10341, 
'Margen': 10341, 
'Municipio': 10341, 
'Precio Biodiesel': 47, 
'Precio Bioetanol': 6, 
'Precio Gas Natural Comprimido': 62, 
'Precio Gas Natural Licuado': 35, 
'Precio Gases licuados del petróleo': 631, 
'Precio Gasoleo A': 10199,   DIESEL
'Precio Gasoleo B': 2385,    OMITIR
'Precio Gasolina 95 Protección': 9866, GASOLINA
'Precio Gasolina  98': 6294,  GASOLINA 98
'Precio Nuevo Gasoleo A': 7266, DIESEL
'Provincia': 10341, 
'Remisión': 10341, 
'Rótulo': 10341,
 'Tipo Venta': 10341, 
 '% BioEtanol': 10341, 
 '% Éster metílico': 10341, 
 'IDEESS': 10341, 'IDMunicipio': 10341, 'IDProvincia': 10341, 'IDCCAA': 10341}
'''