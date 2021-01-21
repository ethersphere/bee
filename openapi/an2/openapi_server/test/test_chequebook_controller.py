# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.cheque_all_peers_response import ChequeAllPeersResponse  # noqa: E501
from openapi_server.models.cheque_peer_response import ChequePeerResponse  # noqa: E501
from openapi_server.models.chequebook_address import ChequebookAddress  # noqa: E501
from openapi_server.models.chequebook_balance import ChequebookBalance  # noqa: E501
from openapi_server.models.swap_cashout_status import SwapCashoutStatus  # noqa: E501
from openapi_server.models.transaction_response import TransactionResponse  # noqa: E501
from openapi_server.test import BaseTestCase


class TestChequebookController(BaseTestCase):
    """ChequebookController integration test stubs"""

    def test_chequebook_address_get(self):
        """Test case for chequebook_address_get

        Get the address of the chequebook contract used
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/address',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_balance_get(self):
        """Test case for chequebook_balance_get

        Get the balance of the chequebook
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/balance',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_cashout_peer_id_get(self):
        """Test case for chequebook_cashout_peer_id_get

        Get last cashout action for the peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/cashout/{peer_id}'.format(peer_id='peer_id_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_cashout_peer_id_post(self):
        """Test case for chequebook_cashout_peer_id_post

        Cashout the last cheque for the peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/cashout/{peer_id}'.format(peer_id='peer_id_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_cheque_get(self):
        """Test case for chequebook_cheque_get

        Get last cheques for all peers
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/cheque',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_cheque_peer_id_get(self):
        """Test case for chequebook_cheque_peer_id_get

        Get last cheques for the peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/cheque/{peer_id}'.format(peer_id='peer_id_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_deposit_post(self):
        """Test case for chequebook_deposit_post

        Deposit tokens from overlay address into chequebook
        """
        query_string = [('amount', 56)]
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/deposit',
            method='POST',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chequebook_withdraw_post(self):
        """Test case for chequebook_withdraw_post

        Withdraw tokens from the chequebook to the overlay address
        """
        query_string = [('amount', 56)]
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chequebook/withdraw',
            method='POST',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
