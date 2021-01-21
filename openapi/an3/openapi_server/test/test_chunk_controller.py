# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.response import Response  # noqa: E501
from openapi_server.test import BaseTestCase


class TestChunkController(BaseTestCase):
    """ChunkController integration test stubs"""

    def test_chunks_address_delete(self):
        """Test case for chunks_address_delete

        Delete a chunk from local storage
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chunks/{address}'.format(address='address_example'),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chunks_address_get(self):
        """Test case for chunks_address_get

        Check if chunk at address exists locally
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/chunks/{address}'.format(address='address_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
