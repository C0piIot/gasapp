from django.conf import settings

def build_version(request):
    return {
        "BUILD_VERSION": settings.BUILD_VERSION
    }