import connexion
import six

from openapi_server.models.address import Address  # noqa: E501
from openapi_server.models.new_tag_request import NewTagRequest  # noqa: E501
from openapi_server.models.new_tag_response import NewTagResponse  # noqa: E501
from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.tags_list import TagsList  # noqa: E501
from openapi_server import util


def tags_get():  # noqa: E501
    """Get list of tags

     # noqa: E501


    :rtype: TagsList
    """
    return 'do some magic!'


def tags_post(new_tag_request):  # noqa: E501
    """Create Tag

     # noqa: E501

    :param new_tag_request: 
    :type new_tag_request: dict | bytes

    :rtype: NewTagResponse
    """
    if connexion.request.is_json:
        new_tag_request = NewTagRequest.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'


def tags_uid_delete(uid):  # noqa: E501
    """Delete Tag information using Uid

     # noqa: E501

    :param uid: Uid
    :type uid: int

    :rtype: None
    """
    return 'do some magic!'


def tags_uid_get(uid):  # noqa: E501
    """Get Tag information using Uid

     # noqa: E501

    :param uid: Uid
    :type uid: int

    :rtype: NewTagResponse
    """
    return 'do some magic!'


def tags_uid_patch(uid, address=None):  # noqa: E501
    """Update Total Count and swarm hash for a tag of an input stream of unknown size using Uid

     # noqa: E501

    :param uid: Uid
    :type uid: int
    :param address: Can contain swarm hash to use for the tag
    :type address: dict | bytes

    :rtype: Status
    """
    if connexion.request.is_json:
        address = Address.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'
