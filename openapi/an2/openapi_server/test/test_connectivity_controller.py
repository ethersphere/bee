# coding: utf-8

from __future__ import absolute_import
import unittest

from flask import json
from six import BytesIO

from openapi_server.models.address import Address  # noqa: E501
from openapi_server.models.addresses import Addresses  # noqa: E501
from openapi_server.models.bzz_topology import BzzTopology  # noqa: E501
from openapi_server.models.peers import Peers  # noqa: E501
from openapi_server.models.response import Response  # noqa: E501
from openapi_server.models.rtt_ms import RttMs  # noqa: E501
from openapi_server.models.status import Status  # noqa: E501
from openapi_server.models.welcome_message import WelcomeMessage  # noqa: E501
from openapi_server.test import BaseTestCase


class TestConnectivityController(BaseTestCase):
    """ConnectivityController integration test stubs"""

    def test_addresses_get(self):
        """Test case for addresses_get

        Get overlay and underlay addresses of the node
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/addresses',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_blocklist_get(self):
        """Test case for blocklist_get

        Get a list of blocklisted peers
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/blocklist',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_connect_multi_address_post(self):
        """Test case for connect_multi_address_post

        Connect to address
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/connect/{multi_address}'.format(multi_address='multi_address_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_peers_address_delete(self):
        """Test case for peers_address_delete

        Remove peer
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/peers/{address}'.format(address='address_example'),
            method='DELETE',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_peers_get(self):
        """Test case for peers_get

        Get a list of peers
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/peers',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_pingpong_peer_id_post(self):
        """Test case for pingpong_peer_id_post

        Try connection to node
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/pingpong/{peer_id}'.format(peer_id='peer_id_example'),
            method='POST',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_topology_get(self):
        """Test case for topology_get

        
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/topology',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_welcome_message_get(self):
        """Test case for welcome_message_get

        Get configured P2P welcome message
        """
        headers = { 
            'Accept': 'application/json',
        }
        response = self.client.open(
            '/welcome-message',
            method='GET',
            headers=headers)
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))

    def test_welcome_message_post(self):
        """Test case for welcome_message_post

        Set P2P welcome message
        """
        welcome_message = {
  "welcome_message" : "welcome_message"
}
        headers = { 
            'Accept': 'application/json',
            'Content-Type': 'application/json',
        }
        response = self.client.open(
            '/welcome-message',
            method='POST',
            headers=headers,
            data=json.dumps(welcome_message),
            content_type='application/json')
        self.assert200(response,
                       'Response body is : ' + response.data.decode('utf-8'))


if __name__ == '__main__':
    unittest.main()
