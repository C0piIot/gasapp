from django.core.management.base import BaseCommand
from stations.models import Station

class Command(BaseCommand):
    help = 'Update stations pricing'
    
    def handle(self, *args, **options):
        Station.update_prices()