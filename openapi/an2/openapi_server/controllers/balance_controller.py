import connexion
import six

from openapi_server.models.balance import Balance  # noqa: E501
from openapi_server.models.balances import Balances  # noqa: E501
from openapi_server import util


def balances_address_get(address):  # noqa: E501
    """Get the balances with a specific peer including prepaid services

     # noqa: E501

    :param address: Swarm address of peer
    :type address: str

    :rtype: Balance
    """
    return 'do some magic!'


def balances_get():  # noqa: E501
    """Get the balances with all known peers including prepaid services

     # noqa: E501


    :rtype: Balances
    """
    return 'do some magic!'


def consumed_address_get(address):  # noqa: E501
    """Get the past due consumption balance with a specific peer

     # noqa: E501

    :param address: Swarm address of peer
    :type address: str

    :rtype: Balance
    """
    return 'do some magic!'


def consumed_get():  # noqa: E501
    """Get the past due consumption balances with all known peers

     # noqa: E501


    :rtype: Balances
    """
    return 'do some magic!'
