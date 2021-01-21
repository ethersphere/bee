# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.new_tag_debug_response import NewTagDebugResponse  # noqa: E501
from openapi_server.test import BaseTestCase


class TestTagController(BaseTestCase):
    """TagController integration test stubs"""

    def test_tags_uid_get(self):
        """Test case for tags_uid_get

        Get Tag information using Uid
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/tags/{uid}'.format(uid=56),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
