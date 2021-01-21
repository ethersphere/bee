# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.test import BaseTestCase


class TestSingleOwnerChunkController(BaseTestCase):
    """SingleOwnerChunkController integration test stubs"""

    def test_soc_owner_id_post(self):
        """Test case for soc_owner_id_post

        Upload single owner chunk
        """
        query_string = [('sig', 'sig_example')]
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/soc/{owner}/{id}'.format(owner='owner_example', id='id_example'),
            method='POST',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
