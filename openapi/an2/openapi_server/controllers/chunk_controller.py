import connexion
import six

from openapi_server.models.response import Response  # noqa: E501
from openapi_server import util


def chunks_address_delete(address):  # noqa: E501
    """Delete a chunk from local storage

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def chunks_address_get(address):  # noqa: E501
    """Check if chunk at address exists locally

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'
