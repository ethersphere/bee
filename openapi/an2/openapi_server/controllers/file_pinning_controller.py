import connexion
import six

from openapi_server.models.response import Response  # noqa: E501
from openapi_server import util


def pin_files_address_delete(address):  # noqa: E501
    """Unpin file chunks with given address

     # noqa: E501

    :param address: Swarm address of the file
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def pin_files_address_post(address):  # noqa: E501
    """Pin file with given address

     # noqa: E501

    :param address: Swarm address of the file
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'
