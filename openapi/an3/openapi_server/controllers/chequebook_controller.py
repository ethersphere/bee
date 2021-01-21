import connexion
import six

from openapi_server.models.cheque_all_peers_response import ChequeAllPeersResponse  # noqa: E501
from openapi_server.models.cheque_peer_response import ChequePeerResponse  # noqa: E501
from openapi_server.models.chequebook_address import ChequebookAddress  # noqa: E501
from openapi_server.models.chequebook_balance import ChequebookBalance  # noqa: E501
from openapi_server.models.swap_cashout_status import SwapCashoutStatus  # noqa: E501
from openapi_server.models.transaction_response import TransactionResponse  # noqa: E501
from openapi_server import util


def chequebook_address_get():  # noqa: E501
    """Get the address of the chequebook contract used

     # noqa: E501


    :rtype: ChequebookAddress
    """
    return 'do some magic!'


def chequebook_balance_get():  # noqa: E501
    """Get the balance of the chequebook

     # noqa: E501


    :rtype: ChequebookBalance
    """
    return 'do some magic!'


def chequebook_cashout_peer_id_get(peer_id):  # noqa: E501
    """Get last cashout action for the peer

     # noqa: E501

    :param peer_id: Swarm address of peer
    :type peer_id: str

    :rtype: SwapCashoutStatus
    """
    return 'do some magic!'


def chequebook_cashout_peer_id_post(peer_id):  # noqa: E501
    """Cashout the last cheque for the peer

     # noqa: E501

    :param peer_id: Swarm address of peer
    :type peer_id: str

    :rtype: TransactionResponse
    """
    return 'do some magic!'


def chequebook_cheque_get():  # noqa: E501
    """Get last cheques for all peers

     # noqa: E501


    :rtype: ChequeAllPeersResponse
    """
    return 'do some magic!'


def chequebook_cheque_peer_id_get(peer_id):  # noqa: E501
    """Get last cheques for the peer

     # noqa: E501

    :param peer_id: Swarm address of peer
    :type peer_id: str

    :rtype: ChequePeerResponse
    """
    return 'do some magic!'


def chequebook_deposit_post(amount):  # noqa: E501
    """Deposit tokens from overlay address into chequebook

     # noqa: E501

    :param amount: amount of tokens to deposit
    :type amount: int

    :rtype: TransactionResponse
    """
    return 'do some magic!'


def chequebook_withdraw_post(amount):  # noqa: E501
    """Withdraw tokens from the chequebook to the overlay address

     # noqa: E501

    :param amount: amount of tokens to withdraw
    :type amount: int

    :rtype: TransactionResponse
    """
    return 'do some magic!'
