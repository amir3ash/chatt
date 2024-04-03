package ws

type TestAuthz struct{}

// WhoCanReadTopic implements whoCanReadTopic.
func (t *TestAuthz) WhoCanReadTopic(topicId string) ([]string, error) {
	return []string{"343", "673"}, nil
}

func (t *TestAuthz) TopicsWhichUserCanRead(userId string, topics []string) (topicIds []string, err error) {
	return topics, nil
}
