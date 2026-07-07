from tools.docsgen.extras import parse_labels

FIXTURE = """\
=Lumbridge,3239,3233,1
=Kingdom Of/Misthalin,3217,3321,2
=Toll/Gate,3278,3227,0
"""


def test_parse_labels():
    labels = parse_labels(FIXTURE)
    assert labels[0] == ("Lumbridge", 3239, 3233, 1)
    assert labels[1] == ("Kingdom Of Misthalin", 3217, 3321, 2)
    assert len(labels) == 3


def test_parse_labels_ignores_blank_and_malformed():
    assert parse_labels("\nnonsense\n=OnlyName\n") == []
