import connexion
import six

from openapi_server.models.status import Status  # noqa: E501
from openapi_server import util


def health_get():  # noqa: E501
    """Get health of node

     # noqa: E501


    :rtype: Status
    """
    return 'do some magic!'


def readiness_get():  # noqa: E501
    """Get readiness state of node

     # noqa: E501


    :rtype: Status
    """
    return 'do some magic!'
