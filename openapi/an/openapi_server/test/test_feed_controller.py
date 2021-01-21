# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.reference_response import ReferenceResponse  # noqa: E501
from openapi_server.test import BaseTestCase


class TestFeedController(BaseTestCase):
    """FeedController integration test stubs"""

    def test_feed_owner_topic_get(self):
        """Test case for feed_owner_topic_get

        Find feed update
        """
        query_string = [('index', 'index_example'),
                        ('at', 56),
                        ('type', 'type_example')]
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/feed/{owner}/{topic}'.format(owner='owner_example', topic='topic_example'),
            method='GET',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_feed_owner_topic_post(self):
        """Test case for feed_owner_topic_post

        Create an initial feed root manifest
        """
        query_string = [('type', 'type_example')]
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/v1/feed/{owner}/{topic}'.format(owner='owner_example', topic='topic_example'),
            method='POST',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
