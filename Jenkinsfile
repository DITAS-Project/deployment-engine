//Jenkins file for the present dockerfile
//WIP = work in progress
pipeline {
  agent any
  stages {
    stage('Build') {
      steps {
        sh 'echo "Building!"'
      }
    }
    stage('Deploy') {
      steps {
        echo 'Deploying!'
      }
    }
  }
  post {
    always {
      echo 'Post action fired'      
    }    
  }
}

