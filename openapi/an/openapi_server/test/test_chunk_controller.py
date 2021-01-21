# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server.test import BaseTestCase


class TestChunkController(BaseTestCase):
    """ChunkController integration test stubs"""

    @unittest.skip("application/octet-stream not supported by Connexion")
    def test_chunks_post(self):
        """Test case for chunks_post

        Upload Chunk
        """
        body = (BytesIO(b'some file data'), 'file.txt')
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'application/octet-stream',
            'swarm_tag': 56,
            'swarm_pin': True,
        }
        response = self.client.open(
            '/v1/chunks',
            method='POST',
            headers=headers,
            data=json.dumps(body),
            content_type='application/octet-stream')
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_chunks_reference_get(self):
        """Test case for chunks_reference_get

        Get Chunk
        """
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/chunks/{reference}'.format(reference={}),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
