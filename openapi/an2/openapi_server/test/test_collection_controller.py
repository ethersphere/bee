# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server.test import BaseTestCase


class TestCollectionController(BaseTestCase):
    """CollectionController integration test stubs"""

    def test_bzz_reference_get(self):
        """Test case for bzz_reference_get

        Get index document from a collection of files
        """
        query_string = [('targets', 'targets_example')]
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/bzz/{reference}'.format(reference={}),
            method='GET',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_bzz_reference_path_get(self):
        """Test case for bzz_reference_path_get

        Get referenced file from a collection of files
        """
        query_string = [('targets', 'targets_example')]
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/bzz/{reference}/{path}'.format(reference={}, path='path_example'),
            method='GET',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    @unittest.skip("application/x-tar not supported by Connexion")
    def test_dirs_post(self):
        """Test case for dirs_post

        Upload a collection
        """
        body = (BytesIO(b'some file data'), 'file.txt')
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'application/x-tar',
            'swarm_tag': 56,
            'swarm_pin': True,
            'swarm_encrypt': True,
            'swarm_index_document': index.html,
            'swarm_error_document': error.html,
        }
        response = self.client.open(
            '/v1/dirs',
            method='POST',
            headers=headers,
            data=json.dumps(body),
            content_type='application/x-tar')
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
