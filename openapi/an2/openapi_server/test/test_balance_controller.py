# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.balance import Balance  # noqa: E501
from openapi_server.models.balances import Balances  # noqa: E501
from openapi_server.test import BaseTestCase


class TestBalanceController(BaseTestCase):
    """BalanceController integration test stubs"""

    def test_balances_address_get(self):
        """Test case for balances_address_get

        Get the balances with a specific peer including prepaid services
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/balances/{address}'.format(address='address_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_balances_get(self):
        """Test case for balances_get

        Get the balances with all known peers including prepaid services
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/balances',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_consumed_address_get(self):
        """Test case for consumed_address_get

        Get the past due consumption balance with a specific peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/consumed/{address}'.format(address='address_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_consumed_get(self):
        """Test case for consumed_get

        Get the past due consumption balances with all known peers
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/consumed',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
