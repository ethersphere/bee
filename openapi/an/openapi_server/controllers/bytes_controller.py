import connexion
import six

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server import util


def bytes_post(swarm_tag=None, swarm_pin=None, swarm_encrypt=None, body=None):  # noqa: E501
    """Upload data

     # noqa: E501

    :param swarm_tag: Associate upload with an existing Tag UID
    :type swarm_tag: int
    :param swarm_pin: Represents the pinning state of the bytes
    :type swarm_pin: bool
    :param swarm_encrypt: Represents the encrypting state of the bytes
    :type swarm_encrypt: bool
    :param body: 
    :type body: str

    :rtype: ReferenceResponse
    """
    return 'do some magic!'


def bytes_reference_get(reference):  # noqa: E501
    """Get referenced data

     # noqa: E501

    :param reference: Swarm address reference to content
    :type reference: dict | bytes

    :rtype: file
    """
    if connexion.request.is_json:
        reference =  SwarmReference.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'
