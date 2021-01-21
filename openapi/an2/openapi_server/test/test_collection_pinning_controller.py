# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.response import Response  # noqa: E501
from openapi_server.test import BaseTestCase


class TestCollectionPinningController(BaseTestCase):
    """CollectionPinningController integration test stubs"""

    def test_pin_bzz_address_delete(self):
        """Test case for pin_bzz_address_delete

        Unpin file chunks with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/bzz/{address}'.format(address='address_example'),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_bzz_address_post(self):
        """Test case for pin_bzz_address_post

        Pin collection with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/bzz/{address}'.format(address='address_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
