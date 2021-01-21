# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.settlement import Settlement  # noqa: E501
from openapi_server.models.settlements import Settlements  # noqa: E501
from openapi_server.test import BaseTestCase


class TestSettlementsController(BaseTestCase):
    """SettlementsController integration test stubs"""

    def test_settlements_address_get(self):
        """Test case for settlements_address_get

        Get amount of sent and received from settlements with a peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/settlements/{address}'.format(address='address_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_settlements_get(self):
        """Test case for settlements_get

        Get settlements with all known peers and total amount sent or received
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/settlements',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
