# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.models.swarm_reference import SwarmReference  # noqa: E501
from openapi_server.test import BaseTestCase


class TestFileController(BaseTestCase):
    """FileController integration test stubs"""

    @unittest.skip("Connexion does not support multiple consummes. See https://github.com/zalando/connexion/pull/760")
    def test_files_post(self):
        """Test case for files_post

        Upload file
        """
        query_string = [('name', 'name_example')]
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'multipart/form-data',
            'swarm_tag': 56,
            'swarm_pin': True,
            'swarm_encrypt': True,
            'content_type': 'content_type_example',
        }
        data = dict(file=(BytesIO(b'some file data'), 'file.txt'))
        response = self.client.open(
            '/v1/files',
            method='POST',
            headers=headers,
            data=data,
            content_type='multipart/form-data',
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_files_reference_get(self):
        """Test case for files_reference_get

        Get referenced file
        """
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/files/{reference}'.format(reference={}),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
