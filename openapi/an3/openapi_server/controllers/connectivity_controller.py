import connexion
import six

from openapi_server.models.address import Address  # noqa: E501
from openapi_server.models.addresses import Addresses  # noqa: E501
from openapi_server.models.bzz_topology import BzzTopology  # noqa: E501
from openapi_server.models.peers import Peers  # noqa: E501
from openapi_server.models.response import Response  # noqa: E501
from openapi_server.models.rtt_ms import RttMs  # noqa: E501
from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.welcome_message import WelcomeMessage  # noqa: E501
from openapi_server import util


def addresses_get():  # noqa: E501
    """Get overlay and underlay addresses of the node

     # noqa: E501


    :rtype: Addresses
    """
    return 'do some magic!'


def blocklist_get():  # noqa: E501
    """Get a list of blocklisted peers

     # noqa: E501


    :rtype: Peers
    """
    return 'do some magic!'


def connect_multi_address_post(multi_address):  # noqa: E501
    """Connect to address

     # noqa: E501

    :param multi_address: Underlay address of peer
    :type multi_address: str

    :rtype: Address
    """
    return 'do some magic!'


def peers_address_delete(address):  # noqa: E501
    """Remove peer

     # noqa: E501

    :param address: Swarm address of peer
    :type address: str

    :rtype: Response
    """
    return 'do some magic!'


def peers_get():  # noqa: E501
    """Get a list of peers

     # noqa: E501


    :rtype: Peers
    """
    return 'do some magic!'


def pingpong_peer_id_post(peer_id):  # noqa: E501
    """Try connection to node

     # noqa: E501

    :param peer_id: Swarm address of peer
    :type peer_id: str

    :rtype: RttMs
    """
    return 'do some magic!'


def topology_get():  # noqa: E501
    """topology_get

    Get topology of known network # noqa: E501


    :rtype: BzzTopology
    """
    return 'do some magic!'


def welcome_message_get():  # noqa: E501
    """Get configured P2P welcome message

     # noqa: E501


    :rtype: WelcomeMessage
    """
    return 'do some magic!'


def welcome_message_post(welcome_message=None):  # noqa: E501
    """Set P2P welcome message

     # noqa: E501

    :param welcome_message: 
    :type welcome_message: dict | bytes

    :rtype: Status
    """
    if connexion.request.is_json:
        welcome_message = WelcomeMessage.from_dict(connexion.request.get_json())  # noqa: E501
    return 'do some magic!'
