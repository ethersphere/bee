import connexion
import six

from openapi_server.models.settlement import Settlement  # noqa: E501
from openapi_server.models.settlements import Settlements  # noqa: E501
from openapi_server import util


def settlements_address_get(address):  # noqa: E501
    """Get amount of sent and received from settlements with a peer

     # noqa: E501

    :param address: Swarm address of peer
    :type address: str

    :rtype: Settlement
    """
    return 'do some magic!'


def settlements_get():  # noqa: E501
    """Get settlements with all known peers and total amount sent or received

     # noqa: E501


    :rtype: Settlements
    """
    return 'do some magic!'
