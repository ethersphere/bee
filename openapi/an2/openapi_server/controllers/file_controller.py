import connexion
import six

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server import util


def files_post(name=None, swarm_tag=None, swarm_pin=None, swarm_encrypt=None, content_type=None, file=None):  # noqa: E501
    """Upload file

     # noqa: E501

    :param name: Filename
    :type name: str
    :param swarm_tag: Associate upload with an existing Tag UID
    :type swarm_tag: int
    :param swarm_pin: Represents the pinning state of the file
    :type swarm_pin: bool
    :param swarm_encrypt: Represents the encrypting state of the file
    :type swarm_encrypt: bool
    :param content_type: 
    :type content_type: str
    :param file: 
    :type file: List[str]

    :rtype: ReferenceResponse
    """
    return 'do some magic!'


def files_reference_get(reference):  # noqa: E501
    """Get referenced file

     # noqa: E501

    :param reference: Swarm address of content
    :type reference: dict | bytes

    :rtype: file
    """
    if connexion.request.is_json:
        reference =  SwarmReference.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'
