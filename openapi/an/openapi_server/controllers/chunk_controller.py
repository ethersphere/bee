import connexion
import six

from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server import util


def chunks_post(swarm_tag=None, swarm_pin=None, body=None):  # noqa: E501
    """Upload Chunk

     # noqa: E501

    :param swarm_tag: Associate upload with an existing Tag UID
    :type swarm_tag: int
    :param swarm_pin: Represents the pinning state of the chunk
    :type swarm_pin: bool
    :param body: 
    :type body: str

    :rtype: Status
    """
    return 'do some magic!'


def chunks_reference_get(reference):  # noqa: E501
    """Get Chunk

     # noqa: E501

    :param reference: Swarm address of chunk
    :type reference: dict | bytes

    :rtype: file
    """
    if connexion.request.is_json:
        reference =  SwarmReference.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'
