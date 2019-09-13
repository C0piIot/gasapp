# Generated by Django 2.2.5 on 2019-09-12 12:11

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('stations', '0002_auto_20190912_0804'),
    ]

    operations = [
        migrations.RemoveField(
            model_name='station',
            name='petrol',
        ),
        migrations.AddField(
            model_name='station',
            name='petrol95',
            field=models.DecimalField(blank=True, decimal_places=3, max_digits=6, null=True, verbose_name='gasolina 95'),
        ),
        migrations.AddField(
            model_name='station',
            name='petrol98',
            field=models.DecimalField(blank=True, decimal_places=3, max_digits=6, null=True, verbose_name='gasolina 98'),
        ),
    ]
