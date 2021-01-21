import connexion
import six

from openapi_server import util


def pss_send_topic_targets_post(topic, targets, recipient=None):  # noqa: E501
    """Send to recipient or target with Postal Service for Swarm

     # noqa: E501

    :param topic: Topic name
    :type topic: str
    :param targets: Target message address prefix. If multiple targets are specified, only one would be matched.
    :type targets: str
    :param recipient: Recipient publickey
    :type recipient: str

    :rtype: None
    """
    return 'do some magic!'
