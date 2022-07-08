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
    glp = models.DecimalField(_('GLP'), max_digits=6, decimal_places=3, blank=True, null=True)
    location = gis_models.PointField(_('location'))

    class Meta:
        verbose_name = _('station')
        verbose_name_plural = _('stations')
