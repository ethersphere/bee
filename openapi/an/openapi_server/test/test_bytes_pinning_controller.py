# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.response import Response  # noqa: E501
from openapi_server.test import BaseTestCase


class TestBytesPinningController(BaseTestCase):
    """BytesPinningController integration test stubs"""

    def test_pin_bytes_address_delete(self):
        """Test case for pin_bytes_address_delete

        Unpin bytes chunks with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/bytes/{address}'.format(address='address_example'),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_bytes_address_post(self):
        """Test case for pin_bytes_address_post

        Pin bytes with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/bytes/{address}'.format(address='address_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
