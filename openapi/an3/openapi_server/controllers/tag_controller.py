import connexion
import six

from openapi_server.models.new_tag_debug_response import NewTagDebugResponse  # noqa: E501
from openapi_server import util


def tags_uid_get(uid):  # noqa: E501
    """Get Tag information using Uid

     # noqa: E501

    :param uid: Uid
    :type uid: int

    :rtype: NewTagDebugResponse
    """
    return 'do some magic!'
