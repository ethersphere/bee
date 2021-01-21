# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.bzz_chunks_pinned import BzzChunksPinned  # noqa: E501
from openapi_server.models.pinning_state import PinningState  # noqa: E501
from openapi_server.models.response import Response  # noqa: E501
from openapi_server.test import BaseTestCase


class TestChunkPinningController(BaseTestCase):
    """ChunkPinningController integration test stubs"""

    def test_pin_chunks_address_delete(self):
        """Test case for pin_chunks_address_delete

        Unpin chunk with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/chunks/{address}'.format(address='address_example'),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_chunks_address_get(self):
        """Test case for pin_chunks_address_get

        Get pinning status of chunk with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/chunks/{address}'.format(address='address_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_chunks_address_post(self):
        """Test case for pin_chunks_address_post

        Pin chunk with given address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/chunks/{address}'.format(address='address_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_chunks_address_put(self):
        """Test case for pin_chunks_address_put

        Update chunk pin counter
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/chunks/{address}'.format(address='address_example'),
            method='PUT',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pin_chunks_get(self):
        """Test case for pin_chunks_get

        Get list of pinned chunks
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/pin/chunks',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
