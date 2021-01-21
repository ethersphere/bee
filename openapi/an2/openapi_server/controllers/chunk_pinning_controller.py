import connexion
import six

from openapi_server.models.bzz_chunks_pinned import BzzChunksPinned  # noqa: E501
from openapi_server.models.pinning_state import PinningState  # noqa: E501
from openapi_server.models.response import Response  # noqa: E501
from openapi_server import util


def pin_chunks_address_delete(address):  # noqa: E501
    """Unpin chunk with given address

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def pin_chunks_address_get(address):  # noqa: E501
    """Get pinning status of chunk with given address

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: PinningState
    """
    return 'do some magic!'


def pin_chunks_address_post(address):  # noqa: E501
    """Pin chunk with given address

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def pin_chunks_address_put(address):  # noqa: E501
    """Update chunk pin counter

     # noqa: E501

    :param address: Swarm address of chunk
    :type address: str

    :rtype: PinningState
    """
    return 'do some magic!'


def pin_chunks_get():  # noqa: E501
    """Get list of pinned chunks

     # noqa: E501


    :rtype: BzzChunksPinned
    """
    return 'do some magic!'
