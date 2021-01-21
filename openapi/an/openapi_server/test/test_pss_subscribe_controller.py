# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.test import BaseTestCase


class TestPssSubscribeController(BaseTestCase):
    """PssSubscribeController integration test stubs"""

    def test_pss_subscribe_topic_get(self):
        """Test case for pss_subscribe_topic_get

        Subscribe for messages on the given topic.
        """
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/pss/subscribe/{topic}'.format(topic='topic_example'),
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
