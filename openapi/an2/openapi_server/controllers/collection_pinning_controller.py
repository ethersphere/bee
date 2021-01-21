import connexion
import six

from openapi_server.models.response import Response  # noqa: E501
from openapi_server import util


def pin_bzz_address_delete(address):  # noqa: E501
    """Unpin file chunks with given address

     # noqa: E501

    :param address: Swarm address of the collection
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def pin_bzz_address_post(address):  # noqa: E501
    """Pin collection with given address

     # noqa: E501

    :param address: Swarm address of the collection
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'
