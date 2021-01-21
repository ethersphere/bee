# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.address import Address  # noqa: E501
from openapi_server.models.new_tag_request import NewTagRequest  # noqa: E501
from openapi_server.models.new_tag_response import NewTagResponse  # noqa: E501
from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.tags_list import TagsList  # noqa: E501
from openapi_server.test import BaseTestCase


class TestTagController(BaseTestCase):
    """TagController integration test stubs"""

    def test_tags_get(self):
        """Test case for tags_get

        Get list of tags
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/tags',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_tags_post(self):
        """Test case for tags_post

        Create Tag
        """
        new_tag_request = {
  "address" : "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'application/json',
        }
        response = self.client.open(
            '/v1/tags',
            method='POST',
            headers=headers,
            data=json.dumps(new_tag_request),
            content_type='application/json')
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_tags_uid_delete(self):
        """Test case for tags_uid_delete

        Delete Tag information using Uid
        """
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/tags/{uid}'.format(uid=56),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_tags_uid_get(self):
        """Test case for tags_uid_get

        Get Tag information using Uid
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/tags/{uid}'.format(uid=56),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_tags_uid_patch(self):
        """Test case for tags_uid_patch

        Update Total Count and swarm hash for a tag of an input stream of unknown size using Uid
        """
        address = {
  "address" : "36b7efd913ca4cf880b8eeac5093fa27b0825906c600685b6abdd6566e6cfe8f"
}
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'application/json',
        }
        response = self.client.open(
            '/v1/tags/{uid}'.format(uid=56),
            method='PATCH',
            headers=headers,
            data=json.dumps(address),
            content_type='application/json')
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
