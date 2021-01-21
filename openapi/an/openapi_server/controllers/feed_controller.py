import connexion
import six

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server import util


def feeds_owner_topic_get(owner, topic, index=None, at=None, type=None):  # noqa: E501
    """Find feed update

     # noqa: E501

    :param owner: Owner
    :type owner: str
    :param topic: Topic
    :type topic: str
    :param index: Feed update index
    :type index: str
    :param at: Timestamp of the update (default: now)
    :type at: int
    :param type: Feed indexing scheme (default: sequence)
    :type type: str

    :rtype: ReferenceResponse
    """
    return 'do some magic!'


def feeds_owner_topic_post(owner, topic, type=None):  # noqa: E501
    """Create an initial feed root manifest

     # noqa: E501

    :param owner: Owner
    :type owner: str
    :param topic: Topic
    :type topic: str
    :param type: Feed indexing scheme (default: sequence)
    :type type: str

    :rtype: ReferenceResponse
    """
    return 'do some magic!'
