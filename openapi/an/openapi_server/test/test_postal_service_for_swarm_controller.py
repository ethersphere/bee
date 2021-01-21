# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.test import BaseTestCase


class TestPostalServiceForSwarmController(BaseTestCase):
    """PostalServiceForSwarmController integration test stubs"""

    def test_pss_send_topic_targets_post(self):
        """Test case for pss_send_topic_targets_post

        Send to recipient or target with Postal Service for Swarm
        """
        query_string = [('recipient', 'recipient_example')]
        headers = { 
            'Accept': 'application/problem+json',
        }
        response = self.client.open(
            '/v1/pss/send/{topic}/{targets}'.format(topic='topic_example', targets='targets_example'),
            method='POST',
            headers=headers,
            query_string=query_string)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
