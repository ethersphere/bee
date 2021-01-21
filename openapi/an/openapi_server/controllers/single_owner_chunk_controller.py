import connexion
import six

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server import util


def soc_owner_id_post(owner, id, sig):  # noqa: E501
    """Upload single owner chunk

     # noqa: E501

    :param owner: Owner
    :type owner: str
    :param id: Id
    :type id: str
    :param sig: Signature
    :type sig: str

    :rtype: ReferenceResponse
    """
    return 'do some magic!'
