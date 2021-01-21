import connexion
import six

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server import util


def bzz_reference_get(reference, targets=None):  # noqa: E501
    """Get index document from a collection of files

     # noqa: E501

    :param reference: Swarm address of content
    :type reference: dict | bytes
    :param targets: Global pinning targets prefix
    :type targets: str

    :rtype: file
    """
    if connexion.request.is_json:
        reference =  SwarmReference.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'


def bzz_reference_path_get(reference, path, targets=None):  # noqa: E501
    """Get referenced file from a collection of files

     # noqa: E501

    :param reference: Swarm address of content
    :type reference: dict | bytes
    :param path: Path to the file in the collection.
    :type path: str
    :param targets: Global pinning targets prefix
    :type targets: str

    :rtype: file
    """
    if connexion.request.is_json:
        reference =  SwarmReference.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'


def dirs_post(swarm_tag=None, swarm_pin=None, swarm_encrypt=None, swarm_index_document=None, swarm_error_document=None, body=None):  # noqa: E501
    """Upload a collection

     # noqa: E501

    :param swarm_tag: Associate upload with an existing Tag UID
    :type swarm_tag: int
    :param swarm_pin: Represents the pinning state of the collection
    :type swarm_pin: bool
    :param swarm_encrypt: Represents the encrypting state of the collection
    :type swarm_encrypt: bool
    :param swarm_index_document: Default file to be referenced on path, if exists under that path
    :type swarm_index_document: str
    :param swarm_error_document: Configure custom error document to be returned when a specified path can not be found in collection
    :type swarm_error_document: str
    :param body: 
    :type body: str

    :rtype: ReferenceResponse
    """
    return 'do some magic!'
